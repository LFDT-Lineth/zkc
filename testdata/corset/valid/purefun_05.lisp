(defpurefun (eq (x :binary) (y :binary)) (* (- x y) (- x y)))
;;
(defcolumns (X :binary) (Y :binary))
;; X == 1 || X == Y
(defconstraint c1 () (== 0 (* (- X 1) (eq X Y))))
