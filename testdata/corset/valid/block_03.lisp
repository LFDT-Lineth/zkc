(defcolumns (X :i16) (Y :i16) (Z :i16))
(defconstraint c1 ()
  (if (== X 0)
      ;; if X==0 then Y == Z
      (begin
       (== 0 Y)
       (== 0 Z))
      ;; else X == Y && (Y == 0 || Z == 0)
      (begin
       (== 0 (- X Y))
       (== 0 (* Y Z)))))
