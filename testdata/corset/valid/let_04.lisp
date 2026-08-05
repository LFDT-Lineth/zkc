(defcolumns (X :i16) (Y :i16) (Z :i16))
(defconstraint c1 ()
  (let ((XeqY (- X Y)))
    (if (== 0 X)
        ;; if X==0 then Y == Z
        (begin
         (== 0 Y)
         (== 0 Z))
        ;; else X == Y && (Y == 0 || Z == 0)
        (begin
         (== 0 XeqY)
         (== 0 (* Y Z))))))
