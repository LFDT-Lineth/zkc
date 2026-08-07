;;error:6:40-41:expected u1, found u16
;;error:7:40-41:expected u1, found u16
(defpurefun (eq (x :binary) (y :binary)) (* (- x y) (- x y)))
;;
(defcolumns (X :binary) (Y :i16) (Z :i16))
(defconstraint c1 () (== (- X 1) (eq X Y)))
(defconstraint c2 () (== (- X 1) (eq X Z)))
