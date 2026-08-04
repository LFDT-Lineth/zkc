(defcolumns (X :i32 :padding 1) (Y :i32 :padding 1))
;;
(defconstraint c1 ()
  (if (== X 0)
      (== 1 0)
      (== X Y)))
